#!/usr/bin/env -S jennifer run
# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

/**
 * The mikrotik module (modules/mikrotik.j): a RouterOS API client. Log in to a
 * router and print its interfaces and IP addresses. Needs the default `jennifer`
 * binary (net) and a reachable router with the API service enabled. Pass host /
 * user / password as the first three arguments (default 192.168.88.1 /
 * admin / empty) - over the plain API port, so only on a link you trust.
 * Without a router the connect throws, which this demo catches.
 * Run: jennifer run examples/modules/mikrotik_demo.j [host] [user] [password]
 * @module mikrotik_demo
 */
use io;
use os;
import "../../modules/mikrotik.j" as mikrotik;

def host as string init "192.168.88.1";
def user as string init "admin";
def password as string init "";
if (len(os.ARGS) > 1) {
    $host = os.ARGS[1];
}
if (len(os.ARGS) > 2) {
    $user = os.ARGS[2];
}
if (len(os.ARGS) > 3) {
    $password = os.ARGS[3];
}

io.printf("connecting to %s (user %s) ...\n", $host, $user);
try {
    # The plain API port (8728) sends the password in the clear, so connect
    # refuses it unless allowInsecureAuth is set: fine on a lab link, otherwise
    # use mikrotik.optionsTLS (api-ssl, 8729).
    def o as mikrotik.Options init mikrotik.options($host, $user, $password);
    $o.allowInsecureAuth = true;
    def s as mikrotik.Session init mikrotik.connect($o);
    io.printf("logged in\n\ninterfaces:\n");
    for (def iface in mikrotik.print($s, "/interface")) {
        io.printf(
            "  %s|pad=14 %s|pad=10 running=%s\n",
            $iface["name"],
            $iface["type"],
            $iface["running"]);
    }
    io.printf("\nip addresses:\n");
    for (def addr in mikrotik.print($s, "/ip/address")) {
        io.printf("  %s|pad=20 on %s\n", $addr["address"], $addr["interface"]);
    }
    mikrotik.close($s);
} catch (e) {
    io.printf("router unavailable: %s\n", $e.message);
}
