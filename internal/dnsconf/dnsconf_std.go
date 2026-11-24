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
	"strings"
	"sync"
	"time"
)

// The default binary's resolver (M29.5). Go's pure resolver reads only
// /etc/resolv.conf and /etc/hosts and offers no way to name other files, but
// it does take a Dial function. When the chosen files are not the system's,
// Setup sets net.DefaultResolver to PreferGo with a Dial that returns one end
// of an in-process pipe: a stub on the other end reads Go's query, answers it
// from the chosen hosts file (A, AAAA and PTR of a listed name) or forwards
// it byte for byte to the chosen resolv.conf's nameservers, one after the
// other. Every lookup in the process goes through net.DefaultResolver (the
// net library, sql's drivers, every dialer without a resolver of its own), so
// the files apply everywhere.
//
// Go still reads /etc/resolv.conf for the search list, ndots, timeout and
// attempts (its defaults when the file is missing), and /etc/hosts before
// asking the stub; only the nameservers and the hosts entries come from the
// chosen files.

// CustomResolver is true on the default binary, which routes lookups through
// the chosen files; jennifer-tiny has no network library.
const CustomResolver = true

var (
	setupOnce sync.Once
	active    Config
)

// Setup picks the files from the environment once per process and, when they
// are not the system's, routes net.DefaultResolver through the stub. It
// returns the configuration in effect. Safe to call from every interpreter.
func Setup() Config {
	setupOnce.Do(func() {
		active = Load()
		if active.Custom() {
			Install(net.DefaultResolver, active)
		}
	})
	return active
}

// SetForTest makes Setup report c (the process resolver is left as it is) and
// returns the function restoring the previous configuration. For the tests of
// the libraries that explain a failed lookup.
func SetForTest(c Config) func() {
	Setup()
	old := active
	active = c
	return func() { active = old }
}

// Install points r at a stub serving c. Exposed for tests, which use their own
// Resolver instead of the process-wide one.
func Install(r *net.Resolver, c Config) {
	s := &stub{conf: c}
	r.PreferGo = true
	r.Dial = s.dial
}

// stub answers one Go resolver exchange per dial.
type stub struct {
	conf Config
}

// attemptTimeout bounds one exchange with one server when the caller set no
// deadline; resolv.conf's own default `timeout:5`.
const attemptTimeout = 5 * time.Second

func (s *stub) dial(ctx context.Context, network, _ string) (net.Conn, error) {
	if h := s.conf.Hint(); h != "" && (s.conf.Resolv.Missing || s.conf.Hosts.Missing) {
		return nil, errors.New(h)
	}
	servers, hosts, err := s.load()
	if err != nil {
		return nil, err
	}
	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Now().Add(attemptTimeout)
	}
	client, server := net.Pipe()
	tcp := strings.HasPrefix(network, "tcp")
	go serve(server, servers, hosts, tcp, deadline)
	return client, nil
}

// load reads the chosen files on every exchange, so an edit applies to the
// next lookup (Go re-reads /etc/resolv.conf every few seconds too).
func (s *stub) load() ([]string, *hostsTable, error) {
	servers := defaultServers
	if p := s.conf.Resolv.Path; p != "" {
		data, err := os.ReadFile(p)
		if err != nil {
			return nil, nil, err
		}
		if ns := parseResolvConf(data); len(ns) > 0 {
			servers = ns
		}
	}
	var hosts *hostsTable
	if f := s.conf.Hosts; f.Path != "" && (f.Source == SourceEnv || f.Source == SourceBuild) {
		data, err := os.ReadFile(f.Path)
		if err != nil {
			return nil, nil, err
		}
		hosts = parseHosts(data)
	}
	return servers, hosts, nil
}

// serve runs the stub's end of the pipe: Go's resolver speaks the stream form
// (a two-byte length before each message) to a conn that is not a PacketConn.
func serve(c net.Conn, servers []string, hosts *hostsTable, tcp bool, deadline time.Time) {
	defer c.Close()
	_ = c.SetDeadline(deadline)
	for {
		var lb [2]byte
		if _, err := io.ReadFull(c, lb[:]); err != nil {
			return
		}
		query := make([]byte, binary.BigEndian.Uint16(lb[:]))
		if _, err := io.ReadFull(c, query); err != nil {
			return
		}
		resp := answerQuery(query, servers, hosts, tcp, deadline)
		if resp == nil {
			// Every server timed out: hold the pipe open so the client's own
			// deadline fires and Go reports a timeout, not a closed stream.
			time.Sleep(time.Until(deadline))
			return
		}
		out := binary.BigEndian.AppendUint16(make([]byte, 0, 2+len(resp)), uint16(len(resp)))
		if _, err := c.Write(append(out, resp...)); err != nil {
			return
		}
	}
}

// answerQuery answers from the hosts table when it lists the name, else
// forwards. A query that does not parse is forwarded too: the server decides.
func answerQuery(query []byte, servers []string, hosts *hostsTable, tcp bool, deadline time.Time) []byte {
	q, err := parseQuestion(query)
	if err == nil && hosts != nil && q.qclass == classIN {
		if resp := fromHosts(query, q, hosts); resp != nil {
			return resp
		}
	}
	resp, timedOut := forward(query, servers, tcp, deadline)
	if resp == nil && !timedOut && err == nil {
		return reply(query, q, rcodeServFail, 0, nil)
	}
	return resp
}

// fromHosts answers an A / AAAA / PTR question about a name the hosts file
// lists: its addresses of that family (none is an empty, successful answer, as
// for a name the file knows only by its other family), or its names.
func fromHosts(query []byte, q question, hosts *hostsTable) []byte {
	switch q.qtype {
	case typeA, typeAAAA:
		addrs, ok := hosts.byName[q.name]
		if !ok {
			return nil
		}
		var rd [][]byte
		for _, a := range addrs {
			if q.qtype == typeA && a.Is4() {
				b := a.As4()
				rd = append(rd, b[:])
			} else if q.qtype == typeAAAA && a.Is6() {
				b := a.As16()
				rd = append(rd, b[:])
			}
		}
		return reply(query, q, rcodeOK, q.qtype, rd)
	case typePTR:
		names, ok := hosts.byAddr[q.name]
		if !ok {
			return nil
		}
		var rd [][]byte
		for _, n := range names {
			rd = append(rd, appendName(nil, n))
		}
		return reply(query, q, rcodeOK, typePTR, rd)
	}
	return nil
}

// forward relays a query to each server in turn until one answers with
// anything but SERVFAIL / REFUSED, splitting the time left between the
// servers not yet tried. It reports whether the last failure was a timeout.
func forward(query []byte, servers []string, tcp bool, deadline time.Time) ([]byte, bool) {
	var fallback []byte
	timedOut := false
	for i, srv := range servers {
		left := time.Until(deadline)
		if left <= 0 {
			timedOut = true
			break
		}
		until := time.Now().Add(left / time.Duration(len(servers)-i))
		resp, err := exchange(srv, query, tcp, until)
		if err != nil {
			var ne net.Error
			timedOut = errors.As(err, &ne) && ne.Timeout()
			continue
		}
		timedOut = false
		if len(resp) >= 4 {
			if rc := resp[3] & 0xF; rc == rcodeServFail || rc == 5 /* REFUSED */ {
				fallback = resp
				continue
			}
		}
		return resp, false
	}
	if fallback != nil {
		return fallback, false
	}
	return nil, timedOut
}

// exchange sends one message to one server over UDP or TCP and returns the
// reply carrying the same id.
func exchange(server string, msg []byte, tcp bool, deadline time.Time) ([]byte, error) {
	network := "udp"
	if tcp {
		network = "tcp"
	}
	ctx, cancel := context.WithDeadline(context.Background(), deadline)
	defer cancel()
	var d net.Dialer
	c, err := d.DialContext(ctx, network, server)
	if err != nil {
		return nil, err
	}
	defer c.Close()
	_ = c.SetDeadline(deadline)
	id := binary.BigEndian.Uint16(msg)
	if tcp {
		out := binary.BigEndian.AppendUint16(make([]byte, 0, 2+len(msg)), uint16(len(msg)))
		if _, err := c.Write(append(out, msg...)); err != nil {
			return nil, err
		}
		var lb [2]byte
		if _, err := io.ReadFull(c, lb[:]); err != nil {
			return nil, err
		}
		resp := make([]byte, binary.BigEndian.Uint16(lb[:]))
		if _, err := io.ReadFull(c, resp); err != nil {
			return nil, err
		}
		return resp, nil
	}
	if _, err := c.Write(msg); err != nil {
		return nil, err
	}
	buf := make([]byte, 65535)
	for {
		n, err := c.Read(buf)
		if err != nil {
			return nil, err
		}
		if n >= headerLen && binary.BigEndian.Uint16(buf) == id {
			return append([]byte(nil), buf[:n]...), nil
		}
	}
}

// Query looks host up against the named servers only ("ip" or "ip:port"),
// for net.lookupWith's dnsServers: no hosts file, no search list, the name taken as
// absolute. The result is the IPv4 addresses, then the IPv6 ones, in the
// servers' order. A failure is a *net.DNSError like the system resolver's.
func Query(ctx context.Context, servers []string, host string) ([]string, error) {
	if a, err := netip.ParseAddr(host); err == nil {
		return []string{a.String()}, nil
	}
	name := strings.TrimSuffix(host, ".")
	if name == "" || len(name) > 253 {
		return nil, &net.DNSError{Err: "no such host", Name: host, IsNotFound: true}
	}
	for _, label := range strings.Split(name, ".") {
		if label == "" || len(label) > 63 {
			return nil, &net.DNSError{Err: "no such host", Name: host, IsNotFound: true}
		}
	}
	var out []string
	notFound := 0
	var lastErr *net.DNSError
	for i, qtype := range []uint16{typeA, typeAAAA} {
		addrs, err := queryType(ctx, servers, name, qtype, uint16(time.Now().UnixNano())+uint16(i))
		if err != nil {
			if err.IsNotFound {
				notFound++
			}
			lastErr = err
			continue
		}
		for _, a := range addrs {
			out = append(out, a.String())
		}
	}
	if len(out) > 0 {
		return out, nil
	}
	if notFound == 2 || lastErr == nil {
		return nil, &net.DNSError{Err: "no such host", Name: host, IsNotFound: true}
	}
	lastErr.Name = host
	return nil, lastErr
}

// queryType asks each server in turn for one record type: UDP, then TCP when
// the reply is truncated. An NXDOMAIN or an empty answer is IsNotFound.
func queryType(ctx context.Context, servers []string, name string, qtype, id uint16) ([]netip.Addr, *net.DNSError) {
	msg := buildQuery(id, name, qtype)
	var last *net.DNSError
	for _, srv := range servers {
		deadline := time.Now().Add(attemptTimeout)
		if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
			deadline = d
		}
		if ctx.Err() != nil {
			return nil, &net.DNSError{Err: "i/o timeout", Server: srv, IsTimeout: true}
		}
		resp, err := exchange(srv, msg, false, deadline)
		var a answer
		if err == nil {
			a, err = parseReply(resp, id, name, qtype)
			if err == nil && a.truncated {
				if resp, err = exchange(srv, msg, true, deadline); err == nil {
					a, err = parseReply(resp, id, name, qtype)
				}
			}
		}
		if err != nil {
			var ne net.Error
			last = &net.DNSError{Err: err.Error(), Server: srv, IsTimeout: errors.As(err, &ne) && ne.Timeout()}
			continue
		}
		switch a.rcode {
		case rcodeOK:
			if len(a.addrs) == 0 {
				return nil, &net.DNSError{Err: "no such host", Server: srv, IsNotFound: true}
			}
			return a.addrs, nil
		case rcodeNXDomain:
			return nil, &net.DNSError{Err: "no such host", Server: srv, IsNotFound: true}
		default:
			last = &net.DNSError{Err: "server misbehaving", Server: srv, IsTemporary: true}
		}
	}
	if last == nil {
		last = &net.DNSError{Err: "no DNS servers", IsTemporary: true}
	}
	return nil, last
}

// ServerAddr normalises a lookupWith dnsServers entry: an IP address, optionally with a
// port ("192.0.2.1", "192.0.2.1:5353", "::1", "[::1]:5353"); port 53 by
// default. A host name is refused: looking it up would need the resolver.
func ServerAddr(s string) (string, error) {
	if a, err := netip.ParseAddr(s); err == nil {
		return net.JoinHostPort(a.String(), "53"), nil
	}
	ap, err := netip.ParseAddrPort(s)
	if err != nil || ap.Port() == 0 {
		return "", errors.New("a DNS server is an IP address with an optional port (\"192.0.2.1\", \"[2001:db8::1]:5353\"), got \"" + s + "\"")
	}
	return ap.String(), nil
}
