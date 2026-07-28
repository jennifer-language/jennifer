#!/usr/bin/env -S jennifer run
# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

/**
 * The ipnet module (modules/ipnet.j): parse IPv4 / IPv6 addresses and CIDR
 * blocks, render them canonically, and run subnet / allow-list math.
 * Run: jennifer run examples/modules/ipnet_demo.j
 * @module ipnet_demo
 */
use io;
use strings;
import "../../modules/ipnet.j" as ipnet;

# scopeLabel names an address's scope by matching the total Scope enum the
# ipnet module returns (a payload-less variant match over a module enum).
func scopeLabel(a as ipnet.Address) {
    match (ipnet.scope($a)) {
        when Global { return "global"; }
        when Private { return "private"; }
        when Loopback { return "loopback"; }
        when LinkLocal { return "link-local"; }
        when Multicast { return "multicast"; }
        when Unspecified { return "unspecified"; }
        when Reserved { return "reserved"; }
    }
    return "?";
}

# Canonical formatting (RFC 5952 for IPv6).
io.printf("=== canonical addresses ===\n");
for (def s in [
    "192.168.1.1",
    "2001:0db8:0000:0000:0000:0000:0000:0001",
    "::1",
    "fe80::1ff:fe23:4567:890a"
]) {
    io.printf("  %s -> %s\n", $s, ipnet.toString(ipnet.parseAddress($s)));
}

# Subnet facts.
io.printf("=== subnet facts ===\n");
for (def cidr in ["192.168.1.0/24", "203.0.113.128/26", "2001:db8::/32"]) {
    def net as ipnet.Network init ipnet.parse($cidr);
    io.printf(
        "  %s  netmask=%s  broadcast=%s\n",
        ipnet.networkString($net),
        ipnet.toString(ipnet.netmask($net)),
        ipnet.toString(ipnet.broadcast($net)));
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

# Subnetting: split a /24 into /26s, then aggregate the halves back.
io.printf("=== split / aggregate ===\n");
def block as ipnet.Network init ipnet.parse("192.168.1.0/24");
io.printf("  %s has %d addresses; usable %s .. %s\n",
    ipnet.networkString($block),
    ipnet.hostCount($block),
    ipnet.toString(ipnet.firstUsable($block)),
    ipnet.toString(ipnet.lastUsable($block)));
def subs as list of ipnet.Network init ipnet.split($block, 26);
def parts as list of string init [];
for (def s in $subs) {
    $parts[] = ipnet.networkString($s);
}
io.printf("  split /24 -> /26: %s\n", strings.join($parts, ", "));
def merged as list of ipnet.Network init ipnet.aggregate([
    ipnet.parse("10.0.0.0/9"),
    ipnet.parse("10.128.0.0/9")
]);
io.printf("  aggregate 10.0.0.0/9 + 10.128.0.0/9 -> %s\n", ipnet.networkString($merged[0]));

# Classification: one match over the total Scope enum.
io.printf("=== scope ===\n");
for (def a in ["8.8.8.8", "10.0.0.1", "127.0.0.1", "169.254.1.1", "192.0.2.5", "fe80::1", "::1"]) {
    io.printf("  %s -> %s\n", $a, scopeLabel(ipnet.parseAddress($a)));
}
