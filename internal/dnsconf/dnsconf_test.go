// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

//go:build !tinygo

package dnsconf

import (
	"context"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"jennifer-lang.dev/jennifer/internal/version"
)

func withPrefix(t *testing.T, p string) {
	t.Helper()
	old := version.Prefix
	version.Prefix = p
	t.Cleanup(func() { version.Prefix = old })
}

func envOf(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func filesOf(paths ...string) func(string) bool {
	return func(p string) bool {
		for _, x := range paths {
			if x == p {
				return true
			}
		}
		return false
	}
}

func TestChoosePrecedence(t *testing.T) {
	withPrefix(t, "/opt/j")
	all := filesOf("/env/resolv.conf", "/opt/j/etc/resolv.conf", "/etc/resolv.conf", "/opt/j/etc/hosts", "/etc/hosts")

	c := Resolve(envOf(map[string]string{ResolvConfEnv: "/env/resolv.conf"}), all)
	if c.Resolv.Path != "/env/resolv.conf" || c.Resolv.Source != SourceEnv || c.Resolv.Missing {
		t.Errorf("env layer: %+v", c.Resolv)
	}
	if c.Hosts.Path != "/opt/j/etc/hosts" || c.Hosts.Source != SourceBuild {
		t.Errorf("hosts build layer: %+v", c.Hosts)
	}
	if !c.Custom() {
		t.Error("an env / build file is custom")
	}

	c = Resolve(envOf(nil), filesOf("/etc/resolv.conf", "/etc/hosts"))
	if c.Resolv.Path != "/etc/resolv.conf" || c.Resolv.Source != SourceSystem {
		t.Errorf("system layer: %+v", c.Resolv)
	}
	if want := []string{"/opt/j/etc/resolv.conf", "/etc/resolv.conf"}; !reflect.DeepEqual(c.Resolv.Tried, want) {
		t.Errorf("tried %v, want %v", c.Resolv.Tried, want)
	}
	if c.Custom() || c.Hint() != "" {
		t.Errorf("system files: custom %v, hint %q", c.Custom(), c.Hint())
	}

	// An env file that does not exist is reported, never skipped.
	c = Resolve(envOf(map[string]string{HostsEnv: "/missing/hosts"}), all)
	if !c.Hosts.Missing || c.Hosts.Path != "/missing/hosts" || !c.Custom() {
		t.Errorf("missing env file: %+v", c.Hosts)
	}
	if h := c.Hint(); !strings.Contains(h, "JENNIFER_HOSTS names \"/missing/hosts\"") {
		t.Errorf("hint %q", h)
	}
}

// The FHS prefix keeps its configuration in /etc: no second layer, so the
// default build looks at exactly the files Go reads itself.
func TestPrefixUsrIsSystem(t *testing.T) {
	for _, p := range []string{"/usr", "/usr/", "/"} {
		withPrefix(t, p)
		c := Resolve(envOf(nil), filesOf("/etc/resolv.conf"))
		if !reflect.DeepEqual(c.Resolv.Tried, []string{"/etc/resolv.conf"}) || c.Custom() {
			t.Errorf("prefix %q: %+v", p, c.Resolv)
		}
	}
	withPrefix(t, "/opt/jennifer/")
	if got := version.SysconfDir(); got != "/opt/jennifer/etc" {
		t.Errorf("SysconfDir = %q", got)
	}
}

// No resolv.conf anywhere: Go's loopback default stays, and the hint names
// every file looked for and the variable.
func TestHintNoConfig(t *testing.T) {
	withPrefix(t, "/opt/j")
	c := Resolve(envOf(nil), filesOf())
	if c.Resolv.Source != SourceNone || c.Custom() {
		t.Fatalf("no config: %+v", c.Resolv)
	}
	h := c.Hint()
	for _, want := range []string{"JENNIFER_RESOLV_CONF", "/opt/j/etc/resolv.conf", "/etc/resolv.conf", "127.0.0.1:53"} {
		if !strings.Contains(h, want) {
			t.Errorf("hint %q lacks %q", h, want)
		}
	}
	msg := c.Explain("net.lookup: x: no such host")
	if c.Explain(msg) != msg {
		t.Error("Explain appends the hint once")
	}
}

func TestParseFiles(t *testing.T) {
	ns := parseResolvConf([]byte("# c\nsearch lan\nnameserver 192.0.2.1\nnameserver  2001:db8::1 # x\nnameserver bogus\nnameserver\n"))
	if want := []string{"192.0.2.1:53", "[2001:db8::1]:53"}; !reflect.DeepEqual(ns, want) {
		t.Errorf("nameservers %v, want %v", ns, want)
	}
	h := parseHosts([]byte("127.0.0.1 localhost\n10.1.2.3 MyHost.test alias.test. # c\nbad line\nfe80::1%lo x.test\n2001:db8::5 myhost.test\n"))
	if got := h.byName["myhost.test"]; len(got) != 2 || got[0].String() != "10.1.2.3" || got[1].String() != "2001:db8::5" {
		t.Errorf("myhost.test -> %v", got)
	}
	if _, ok := h.byName["x.test"]; ok {
		t.Error("a zoned address is skipped")
	}
	if got := h.byAddr["3.2.1.10.in-addr.arpa"]; !reflect.DeepEqual(got, []string{"myhost.test", "alias.test"}) {
		t.Errorf("PTR -> %v", got)
	}
	if got := reverseName(netip.MustParseAddr("2001:db8::5")); !strings.HasPrefix(got, "5.0.0.0.") || !strings.HasSuffix(got, ".8.b.d.0.1.0.0.2.ip6.arpa") {
		t.Errorf("ip6 reverse %q", got)
	}
}

func TestServerAddr(t *testing.T) {
	for in, want := range map[string]string{
		"192.0.2.1": "192.0.2.1:53", "192.0.2.1:5353": "192.0.2.1:5353",
		"::1": "[::1]:53", "[2001:db8::1]:5353": "[2001:db8::1]:5353",
	} {
		if got, err := ServerAddr(in); err != nil || got != want {
			t.Errorf("ServerAddr(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, bad := range []string{"", "dns.example", "dns.example:53", "192.0.2.1:0", "192.0.2.1:x"} {
		if _, err := ServerAddr(bad); err == nil {
			t.Errorf("ServerAddr(%q) accepted", bad)
		}
	}
}

// fakeDNS is a loopback DNS server on one port, UDP and TCP:
// example.test A 192.0.2.7 / AAAA 2001:db8::7; big.test truncated over UDP,
// A 192.0.2.8 over TCP; fail.test SERVFAIL; anything else NXDOMAIN.
type fakeDNS struct {
	addr    string
	port    string
	mu      sync.Mutex
	queries []string
}

func startFakeDNS(t *testing.T) *fakeDNS {
	t.Helper()
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := pc.LocalAddr().String()
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		pc.Close()
		t.Skipf("no TCP listener on the UDP port: %v", err)
	}
	t.Cleanup(func() { pc.Close(); ln.Close() })
	_, port, _ := net.SplitHostPort(addr)
	f := &fakeDNS{addr: addr, port: port}
	go func() {
		buf := make([]byte, 4096)
		for {
			n, peer, err := pc.ReadFrom(buf)
			if err != nil {
				return
			}
			if resp := f.answer(buf[:n], false); resp != nil {
				_, _ = pc.WriteTo(resp, peer)
			}
		}
	}()
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer c.Close()
				var lb [2]byte
				if _, err := io.ReadFull(c, lb[:]); err != nil {
					return
				}
				q := make([]byte, binary.BigEndian.Uint16(lb[:]))
				if _, err := io.ReadFull(c, q); err != nil {
					return
				}
				resp := f.answer(q, true)
				out := binary.BigEndian.AppendUint16(nil, uint16(len(resp)))
				_, _ = c.Write(append(out, resp...))
			}()
		}
	}()
	return f
}

func (f *fakeDNS) answer(query []byte, tcp bool) []byte {
	q, err := parseQuestion(query)
	if err != nil {
		return nil
	}
	f.mu.Lock()
	f.queries = append(f.queries, q.name)
	f.mu.Unlock()
	ip := func(s string) []byte { return netip.MustParseAddr(s).AsSlice() }
	switch {
	case q.name == "example.test" && q.qtype == typeA:
		return reply(query, q, rcodeOK, typeA, [][]byte{ip("192.0.2.7")})
	case q.name == "example.test" && q.qtype == typeAAAA:
		return reply(query, q, rcodeOK, typeAAAA, [][]byte{ip("2001:db8::7")})
	case q.name == "example.test", q.name == "big.test" && q.qtype != typeA:
		return reply(query, q, rcodeOK, q.qtype, nil)
	case q.name == "big.test" && !tcp:
		r := reply(query, q, rcodeOK, typeA, nil)
		r[2] |= 0x02 // TC
		return r
	case q.name == "big.test":
		return reply(query, q, rcodeOK, typeA, [][]byte{ip("192.0.2.8")})
	case q.name == "fail.test":
		return reply(query, q, rcodeServFail, 0, nil)
	}
	return reply(query, q, rcodeNXDomain, 0, nil)
}

func TestQueryLoopbackFake(t *testing.T) {
	f := startFakeDNS(t)
	ctx := context.Background()
	got, err := Query(ctx, []string{f.addr}, "example.test")
	if err != nil || !reflect.DeepEqual(got, []string{"192.0.2.7", "2001:db8::7"}) {
		t.Errorf("example.test -> %v, %v", got, err)
	}
	if got, err := Query(ctx, []string{f.addr}, "Big.Test."); err != nil || !reflect.DeepEqual(got, []string{"192.0.2.8"}) {
		t.Errorf("big.test (truncated, then TCP) -> %v, %v", got, err)
	}
	var dnsErr *net.DNSError
	if _, err := Query(ctx, []string{f.addr}, "nope.test"); !errors.As(err, &dnsErr) || !dnsErr.IsNotFound {
		t.Errorf("nope.test: %v", err)
	}
	if _, err := Query(ctx, []string{f.addr}, "fail.test"); !errors.As(err, &dnsErr) || dnsErr.IsNotFound || !strings.Contains(err.Error(), "server misbehaving") {
		t.Errorf("fail.test: %v", err)
	}
	if got, err := Query(ctx, nil, "192.0.2.1"); err != nil || !reflect.DeepEqual(got, []string{"192.0.2.1"}) {
		t.Errorf("an IP literal: %v, %v", got, err)
	}
	// A second server is asked when the first does not answer.
	silent, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer silent.Close()
	tctx, cancel := context.WithTimeout(ctx, 300*time.Millisecond)
	defer cancel()
	start := time.Now()
	if _, err := Query(tctx, []string{silent.LocalAddr().String()}, "example.test"); !errors.As(err, &dnsErr) || !dnsErr.IsTimeout {
		t.Errorf("silent server: %v", err)
	}
	if time.Since(start) > 3*time.Second {
		t.Errorf("the deadline was not kept: %v", time.Since(start))
	}
}

// writeFile writes a file in a test dir.
func writeFile(t *testing.T, dir, name, body string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// The stub: Go's own resolver, routed through the chosen resolv.conf (whose
// nameserver is the loopback fake) and hosts file.
func TestStubResolver(t *testing.T) {
	f := startFakeDNS(t)
	old := nameserverPort
	nameserverPort = f.port
	t.Cleanup(func() { nameserverPort = old })

	dir := t.TempDir()
	c := Config{
		Resolv: File{Path: writeFile(t, dir, "resolv.conf", "nameserver 127.0.0.1\n"), Source: SourceEnv, Env: ResolvConfEnv},
		Hosts:  File{Path: writeFile(t, dir, "hosts", "10.1.2.3 jhost.test\n"), Source: SourceEnv, Env: HostsEnv},
	}
	r := &net.Resolver{}
	Install(r, c)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if got, err := r.LookupHost(ctx, "example.test"); err != nil || !containsString(got, "192.0.2.7") {
		t.Errorf("example.test via the stub -> %v, %v", got, err)
	}
	if got, err := r.LookupHost(ctx, "jhost.test"); err != nil || !reflect.DeepEqual(got, []string{"10.1.2.3"}) {
		t.Errorf("jhost.test from the hosts file -> %v, %v", got, err)
	}
	if got, err := r.LookupAddr(ctx, "10.1.2.3"); err != nil || !reflect.DeepEqual(got, []string{"jhost.test."}) {
		t.Errorf("PTR from the hosts file -> %v, %v", got, err)
	}
	if got, err := r.LookupHost(ctx, "big.test"); err != nil || !reflect.DeepEqual(got, []string{"192.0.2.8"}) {
		t.Errorf("big.test (TC, then TCP through the stub) -> %v, %v", got, err)
	}
	var dnsErr *net.DNSError
	if _, err := r.LookupHost(ctx, "nope.test"); !errors.As(err, &dnsErr) || !dnsErr.IsNotFound {
		t.Errorf("nope.test: %v", err)
	}
	f.mu.Lock()
	sawJhost := containsString(f.queries, "jhost.test")
	f.mu.Unlock()
	if sawJhost {
		t.Error("a name the hosts file lists went to the server")
	}
}

// A variable naming a missing file fails every lookup, naming the variable.
func TestStubMissingFile(t *testing.T) {
	c := Config{
		Resolv: File{Path: "/nonexistent/resolv.conf", Source: SourceEnv, Env: ResolvConfEnv, Missing: true},
		Hosts:  File{Source: SourceNone, Env: HostsEnv},
	}
	r := &net.Resolver{}
	Install(r, c)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := r.LookupHost(ctx, "example.test")
	if err == nil || !strings.Contains(err.Error(), "JENNIFER_RESOLV_CONF names \"/nonexistent/resolv.conf\"") {
		t.Errorf("missing resolv.conf: %v", err)
	}
}

// With no explicit file, Setup leaves the process resolver alone.
func TestSystemConfigLeavesDefaultResolver(t *testing.T) {
	c := Config{Resolv: File{Path: "/etc/resolv.conf", Source: SourceSystem}, Hosts: File{Path: "/etc/hosts", Source: SourceSystem}}
	if c.Custom() {
		t.Error("the system files need no stub")
	}
}
