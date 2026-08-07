#!/usr/bin/env -S jennifer run
# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

/**
 * The snmp module (modules/snmp.j) as a server: a self-contained SNMP agent and
 * client in one program. It builds a small MIB, serves it on a loopback UDP
 * socket in a spawned goroutine, then queries itself - no external hardware. A
 * handy simulator for testing an SNMP tool, or a template for exposing your own
 * app's metrics over SNMP.
 * Run: jennifer run examples/modules/snmp_agent_demo.j
 * @module snmp_agent_demo
 */
use io;
use net;
use task;
use channel;
import "../../modules/snmp.j" as snmp;

# The MIB the agent serves: one binding per OID, any SNMP value type.
def a as snmp.Agent init snmp.agent("public", snmp.VERSION2C, [
    snmp.stringVar("1.3.6.1.2.1.1.1.0", "Jennifer SNMP agent"),
    snmp.oidVar("1.3.6.1.2.1.1.2.0", "1.3.6.1.4.1.99999"),
    snmp.varbind("1.3.6.1.2.1.1.3.0", "timeTicks", "", 424242),
    snmp.stringVar("1.3.6.1.2.1.1.5.0", "host-01"),
    snmp.intVar("1.3.6.1.2.1.1.7.0", 72)
]);

# Bind the socket first (so there is no start-up race), then serve on it in a
# spawned goroutine while the main program acts as the client. The stop channel
# shuts the agent down gracefully when we are done.
def sock as net.UDPSocket init net.listenUDP("127.0.0.1:0");
def addr as string init net.address($sock);
def stop as channel of bool init channel.make(1);
def server as task of null init spawn { snmp.serveOn($a, $sock, $stop); };

def c as snmp.Client init snmp.clientWith($addr, "public", snmp.VERSION2C, 2000, 3);

io.printf("GET sysDescr + sysUpTime:\n");
for (def vb in snmp.get($c, ["1.3.6.1.2.1.1.1.0", "1.3.6.1.2.1.1.3.0"])) {
    io.printf("  %s [%s] %s\n", $vb.oid, $vb.type, $vb.value);
}

io.printf("\nWALK the system subtree:\n");
for (def vb in snmp.walk($c, "1.3.6.1.2.1.1")) {
    io.printf("  %s [%s] %s\n", $vb.oid, $vb.type, $vb.value);
}

io.printf("\nSET sysName, then GET it back:\n");
snmp.set($c, [snmp.stringVar("1.3.6.1.2.1.1.5.0", "host-renamed")]);
def after as list of snmp.Varbind init snmp.get($c, ["1.3.6.1.2.1.1.5.0"]);
io.printf("  sysName is now: %s\n", $after[0].value);

# Signal the agent to shut down, then wait for it to finish - a graceful stop.
channel.send($stop, true);
task.wait($server);
net.close($sock);
io.printf("\nagent stopped.\n");
