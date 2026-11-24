// SPDX-License-Identifier: LGPL-3.0-only
// SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

//go:build !tinygo

package netlib_test

import (
	"fmt"
	stdnet "net"
	"strings"
	"testing"

	"jennifer-lang.dev/jennifer/internal/dnsconf"
)

// pickUDPAddr reserves an ephemeral loopback UDP port for a Jennifer listener.
func pickUDPAddr(t *testing.T) string {
	t.Helper()
	p, err := stdnet.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := p.LocalAddr().String()
	_ = p.Close()
	return addr
}

// jenniferDNSFake is a loopback DNS server written in Jennifer: a spawned task
// answers two queries (lookupWith asks A, then AAAA) on a UDP socket,
// example.test A 192.0.2.7 and an empty AAAA answer.
const jenniferDNSFake = `
use io;
use net;
use task;

def sock as net.UDPSocket init net.listenUDP(%q);

func answer(q as bytes) {
    def n as int init len($q);
    def isA as bool init $q[$n - 4] == 0 and $q[$n - 3] == 1;
    def r as bytes;
    $r[] = $q[0];
    $r[] = $q[1];
    $r[] = 129;
    $r[] = 128;
    $r[] = 0;
    $r[] = 1;
    $r[] = 0;
    if ($isA) {
        $r[] = 1;
    } else {
        $r[] = 0;
    }
    for (def i in 0..4) {
        $r[] = 0;
    }
    for (def i in 12..$n) {
        $r[] = $q[$i];
    }
    if ($isA) {
        for (def b in [192, 12, 0, 1, 0, 1, 0, 0, 0, 0, 0, 4, 192, 0, 2, 7]) {
            $r[] = $b;
        }
    }
    return $r;
}

def server as task of int init spawn {
    for (def k in 0..2) {
        def d as net.Datagram init net.recvFrom($sock, 512);
        net.sendTo($sock, $d.peer, answer($d.data));
    }
    return 0;
};

def got as list of string init net.lookupWith("example.test", net.ConnectOptions{dnsServers: [%q], timeoutMs: 5000});
task.wait($server);
net.close($sock);
io.printf("%%v\n", $got);
`

// net.lookupWith with dnsServers asks the named servers only: here a DNS
// fake written in Jennifer on loopback UDP.
func TestLookupWithDNSServersJenniferFake(t *testing.T) {
	addr := pickUDPAddr(t)
	out, err := runProg(t, fmt.Sprintf(jenniferDNSFake, addr, addr))
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if strings.TrimSpace(out) != `["192.0.2.7"]` {
		t.Errorf("lookupWith -> %q", out)
	}
}

// A silent server ends in kind "timeout"; a host name among dnsServers, more
// than 16 of them, or a bad timeoutMs is "runtime" misuse. An IP literal
// answers itself; lookup's result for it equals lookupWith's.
func TestLookupWithDNSServersErrors(t *testing.T) {
	silent, err := stdnet.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer silent.Close()
	out, err := runProg(t, fmt.Sprintf(`
use io;
use net;
try {
    net.lookupWith("example.test", net.ConnectOptions{dnsServers: [%q], timeoutMs: 200});
} catch (e) {
    io.printf("%%s\n", $e.kind);
}
try {
    net.lookupWith("example.test", net.ConnectOptions{dnsServers: ["dns.example"]});
} catch (e) {
    io.printf("%%s: %%s\n", $e.kind, $e.message);
}
try {
    net.lookupWith("example.test", net.ConnectOptions{dnsServers: [%q], timeoutMs: -1});
} catch (e) {
    io.printf("%%s\n", $e.kind);
}
def many as list of string;
for (def i in 0..17) {
    $many[] = "127.0.0.1";
}
try {
    net.lookupWith("example.test", net.ConnectOptions{dnsServers: $many});
} catch (e) {
    io.printf("%%s: %%s\n", $e.kind, $e.message);
}
io.printf("%%v\n", net.lookupWith("192.0.2.1", net.ConnectOptions{dnsServers: ["127.0.0.1"]}));
io.printf("%%v\n", net.lookup("127.0.0.1"));
`, silent.LocalAddr().String(), silent.LocalAddr().String()))
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	want := []string{"timeout", "runtime: net.lookupWith: a DNS server is an IP address", "runtime",
		"runtime: net.lookupWith: net.ConnectOptions.dnsServers names 17 servers, at most 16", `["192.0.2.1"]`, `["127.0.0.1"]`}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != len(want) {
		t.Fatalf("output %q", out)
	}
	for i, w := range want {
		if !strings.HasPrefix(lines[i], w) {
			t.Errorf("line %d = %q, want prefix %q", i, lines[i], w)
		}
	}
}

// With no resolver configuration at all, a failed lookup is a NetError of
// reason "dns" whose message names the files looked for and the variable.
func TestLookupNoConfigHint(t *testing.T) {
	restore := dnsconf.SetForTest(dnsconf.Config{
		Resolv: dnsconf.File{Source: dnsconf.SourceNone, Env: dnsconf.ResolvConfEnv, Tried: []string{"/opt/j/etc/resolv.conf", "/etc/resolv.conf"}},
		Hosts:  dnsconf.File{Source: dnsconf.SourceNone, Env: dnsconf.HostsEnv},
	})
	defer restore()
	out, err := runProg(t, `
use io;
use net;
try {
    net.lookup("nonexistent.invalid");
} catch (e) {
    def ne as net.NetError init $e;
    io.printf("%s %s %s\n", $ne.kind, $ne.reason, $ne.message);
}
`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	for _, want := range []string{"net dns ", "JENNIFER_RESOLV_CONF is not set", "/opt/j/etc/resolv.conf, /etc/resolv.conf"} {
		if !strings.Contains(out, want) {
			t.Errorf("output %q lacks %q", out, want)
		}
	}
}
