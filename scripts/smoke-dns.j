#!/usr/bin/env -S jennifer run
# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>
#
# Name resolution without the network, for the release smoke test of every
# architecture (M29.5, M29.6). A task answers DNS queries on a loopback UDP
# socket (example.test A 192.0.2.7, an empty AAAA answer) and
# `net.lookupWith` with `dnsServers` asks it. With JENNIFER_HOSTS naming a
# file that lists smoke.test, `net.lookup("smoke.test")` also goes through the
# configured resolver's hosts answer. Prints one line per check; any failure throws.

use io;
use net;
use os;
use task;

# reply answers one query: the header and question echoed, plus one A record
# (pointing back at the question's name) when the query asked for A.
func reply(q as bytes) {
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

def sock as net.UDPSocket init net.listenUDP("127.0.0.1:0");
def addr as string init net.address($sock);

def server as task of int init spawn {
    for (def k in 0..2) {
        def d as net.Datagram init net.recvFrom($sock, 512);
        net.sendTo($sock, $d.peer, reply($d.data));
    }
    return 0;
};

def got as list of string init net.lookupWith(
    "example.test",
    net.ConnectOptions{dnsServers: [$addr], timeoutMs: 20000});
task.wait($server);
net.close($sock);
if (len($got) != 1 or $got[0] != "192.0.2.7") {
    throw Error{kind: "smoke", message: "lookupWith did not answer 192.0.2.7"};
}
io.printf("lookupWith: %s\n", $got[0]);

if (os.getEnv("JENNIFER_HOSTS") != "") {
    def hosts as list of string init net.lookup("smoke.test");
    io.printf("hosts: %v\n", $hosts);
}
