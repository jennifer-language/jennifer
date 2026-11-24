// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

//go:build !tinygo

package dnsconf

import (
	"bufio"
	"bytes"
	"net"
	"net/netip"
	"os"
	"strconv"
	"strings"
)

// defaultServers is Go's own choice when a resolv.conf names none or none
// exists: the loopback resolver, never a public one.
var defaultServers = []string{"127.0.0.1:53", "[::1]:53"}

// nameserverPort is the port of a resolv.conf nameserver (the file cannot
// name one); a variable so tests can point it at a loopback fake.
var nameserverPort = "53"

// parseResolvConf returns the `nameserver` addresses of a resolv.conf as
// host:port (port 53). Lines that are not a nameserver with an IP address are
// skipped, as glibc and Go do; search / options lines are Go's concern and
// come from /etc/resolv.conf (see the package doc of the stub).
func parseResolvConf(data []byte) []string {
	var out []string
	sc := bufio.NewScanner(bytes.NewReader(data))
	for sc.Scan() {
		line := sc.Text()
		if i := strings.IndexAny(line, "#;"); i >= 0 {
			line = line[:i]
		}
		f := strings.Fields(line)
		if len(f) < 2 || f[0] != "nameserver" {
			continue
		}
		if addr, err := netip.ParseAddr(f[1]); err == nil {
			out = append(out, net.JoinHostPort(addr.String(), nameserverPort))
		}
	}
	return out
}

// hostsTable is a parsed hosts file: names to addresses, and the reverse
// (in-addr.arpa / ip6.arpa) names to host names.
type hostsTable struct {
	byName map[string][]netip.Addr
	byAddr map[string][]string
	// search is the search list Go's resolver applies before it asks for a
	// name as written (see searchList).
	search []string
}

// parseHosts reads a hosts file: an address, then its names, `#` comments.
// A line whose address does not parse, or that carries an IPv6 zone (which a
// DNS answer cannot express), is skipped, as Go's own reader skips it; so is
// a name that is not a valid DNS name (an empty or over-long label).
func parseHosts(data []byte) *hostsTable {
	t := &hostsTable{byName: map[string][]netip.Addr{}, byAddr: map[string][]string{}}
	sc := bufio.NewScanner(bytes.NewReader(data))
	for sc.Scan() {
		line := sc.Text()
		if i := strings.IndexByte(line, '#'); i >= 0 {
			line = line[:i]
		}
		f := strings.Fields(line)
		if len(f) < 2 {
			continue
		}
		addr, err := netip.ParseAddr(f[0])
		if err != nil || addr.Zone() != "" {
			continue
		}
		addr = addr.Unmap()
		rev := reverseName(addr)
		for _, name := range f[1:] {
			name = strings.ToLower(strings.TrimSuffix(name, "."))
			if !validName(name) {
				continue
			}
			if !containsAddr(t.byName[name], addr) {
				t.byName[name] = append(t.byName[name], addr)
			}
			if !containsString(t.byAddr[rev], name) {
				t.byAddr[rev] = append(t.byAddr[rev], name)
			}
		}
	}
	return t
}

// searchList is the search list Go's resolver reads: the last `search` or
// `domain` line of /etc/resolv.conf, else the domain of the host name. A
// variable so tests can set it.
var searchList = func() []string {
	data, _ := os.ReadFile(systemDir + "/resolv.conf")
	if s := parseSearch(data); s != nil {
		return s
	}
	if hn, err := os.Hostname(); err == nil {
		if i := strings.IndexByte(hn, '.'); i >= 0 && i < len(hn)-1 {
			return []string{strings.ToLower(strings.TrimSuffix(hn[i+1:], "."))}
		}
	}
	return nil
}

// parseSearch returns the domains of a resolv.conf's last `search` or
// `domain` line, lower case without the trailing dot; nil when it has none.
func parseSearch(data []byte) []string {
	var out []string
	sc := bufio.NewScanner(bytes.NewReader(data))
	for sc.Scan() {
		line := sc.Text()
		if i := strings.IndexAny(line, "#;"); i >= 0 {
			line = line[:i]
		}
		f := strings.Fields(line)
		if len(f) < 2 || (f[0] != "search" && f[0] != "domain") {
			continue
		}
		out = out[:0:0]
		for _, d := range f[1:] {
			if d = strings.ToLower(strings.TrimSuffix(d, ".")); d != "" {
				out = append(out, d)
			}
		}
	}
	return out
}

// searchVariant reports whether name is a name the hosts file lists with a
// search domain appended, which Go asks for before the name as written.
func (t *hostsTable) searchVariant(name string) bool {
	for _, d := range t.search {
		if base, ok := strings.CutSuffix(name, "."+d); ok {
			if _, listed := t.byName[base]; listed {
				return true
			}
		}
	}
	return false
}

func containsAddr(list []netip.Addr, a netip.Addr) bool {
	for _, x := range list {
		if x == a {
			return true
		}
	}
	return false
}

func containsString(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// reverseName is the PTR name of an address, lower case, no trailing dot.
func reverseName(a netip.Addr) string {
	const hex = "0123456789abcdef"
	if a.Is4() {
		b := a.As4()
		var sb strings.Builder
		for i := 3; i >= 0; i-- {
			sb.WriteString(strconv.Itoa(int(b[i])))
			sb.WriteByte('.')
		}
		sb.WriteString("in-addr.arpa")
		return sb.String()
	}
	b := a.As16()
	var sb strings.Builder
	for i := 15; i >= 0; i-- {
		sb.WriteByte(hex[b[i]&0xF])
		sb.WriteByte('.')
		sb.WriteByte(hex[b[i]>>4])
		sb.WriteByte('.')
	}
	sb.WriteString("ip6.arpa")
	return sb.String()
}
