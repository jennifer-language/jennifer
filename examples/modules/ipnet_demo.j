#!/usr/bin/env -S jennifer run
# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 mplx <jennifer@mplx.dev>

/**
 * The ipnet module (modules/ipnet.j): parse IPv4 / IPv6 addresses and CIDR
 * blocks, render them canonically, and run subnet / allow-list math.
 * Run: jennifer run examples/modules/ipnet_demo.j
 * @module ipnet_demo
 */
use io;
import "../../modules/ipnet.j" as ipnet;

# Canonical formatting (RFC 5952 for IPv6).
io.printf("=== canonical addresses ===\n");
for (def s in ["192.168.1.1", "2001:0db8:0000:0000:0000:0000:0000:0001", "::1", "fe80::1ff:fe23:4567:890a"]) {
    io.printf("  %s -> %s\n", $s, ipnet.toString(ipnet.parseAddress($s)));
}

# Subnet facts.
io.printf("=== subnet facts ===\n");
for (def cidr in ["192.168.1.0/24", "203.0.113.128/26", "2001:db8::/32"]) {
    def net as ipnet.Network init ipnet.parse($cidr);
    io.printf("  %s  netmask=%s  broadcast=%s\n", ipnet.networkString($net),
        ipnet.toString(ipnet.netmask($net)), ipnet.toString(ipnet.broadcast($net)));
}

# Allow-list check: is a client address inside any allowed CIDR?
io.printf("=== allow-list ===\n");
def allowed as list of ipnet.Network init [];
$allowed[] = ipnet.parse("10.0.0.0/8");
$allowed[] = ipnet.parse("192.168.0.0/16");
$allowed[] = ipnet.parse("2001:db8::/32");

for (def client in ["10.4.5.6", "192.168.1.42", "8.8.8.8", "2001:db8:abcd::1"]) {
    def addr as ipnet.Address init ipnet.parseAddress($client);
    def ok as bool init false;
    for (def net in $allowed) {
        if (ipnet.contains($net, $addr)) {
            $ok = true;
        }
    }
    io.printf("  %s -> %t\n", $client, $ok);
}
