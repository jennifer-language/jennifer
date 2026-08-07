#!/usr/bin/env -S jennifer run
# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

/**
 * The snmp module (modules/snmp.j): an SNMP v1 / v2c manager over UDP. GET a
 * handful of system OIDs and walk the system subtree.
 * Run: jennifer run examples/modules/snmp_demo.j [host] [community] [v1|v2c]
 * (defaults: 127.0.0.1, "public", v2c). Point it at a real agent to see output;
 * against nothing it prints the timeout error and exits.
 * @module snmp_demo
 */
use io;
use os;
import "../../modules/snmp.j" as snmp;

def host as string init "127.0.0.1";
def community as string init "public";
def version as int init snmp.VERSION2C;

if (len(os.ARGS) > 1) {
    $host = os.ARGS[1];
}
if (len(os.ARGS) > 2) {
    $community = os.ARGS[2];
}
if (len(os.ARGS) > 3 and os.ARGS[3] == "v1") {
    $version = snmp.VERSION1;
}

def c as snmp.Client init snmp.clientWith($host + ":161", $community, $version, 3000, 1);

# Standard system-group OIDs (RFC 1213).
def names as map of string to string init {
    "1.3.6.1.2.1.1.1.0": "sysDescr",
    "1.3.6.1.2.1.1.3.0": "sysUpTime",
    "1.3.6.1.2.1.1.5.0": "sysName",
    "1.3.6.1.2.1.1.6.0": "sysLocation"
};

try {
    io.printf("GET system OIDs from %s (%s):\n", $host, $community);
    def keys as list of string init [
        "1.3.6.1.2.1.1.1.0", "1.3.6.1.2.1.1.3.0",
        "1.3.6.1.2.1.1.5.0", "1.3.6.1.2.1.1.6.0"
    ];
    for (def vb in snmp.get($c, $keys)) {
        io.printf("  %s [%s] %s\n", $names[$vb.oid], $vb.type, $vb.value);
    }

    io.printf("\nWALK the system subtree:\n");
    def rows as list of snmp.Varbind init snmp.walk($c, "1.3.6.1.2.1.1");
    io.printf("  %d bindings\n", len($rows));
    for (def vb in $rows) {
        io.printf("  %s [%s] %s\n", $vb.oid, $vb.type, $vb.value);
    }
} catch (e) {
    io.printf("snmp error: %s\n", $e.message);
    io.printf("(point the demo at a reachable agent: snmp_demo.j <host> <community> [v1])\n");
}
