# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 <developer@mplx.eu>
#
# net.j - exercises the M16.2 `net` library. A tiny in-process TCP
# echo: a `spawn`'d server accepts one connection and echoes
# whatever it reads; the main flow connects, sends a message, reads
# it back, and prints it.
#
# Uses `:0` on the server so the kernel picks an ephemeral port,
# then `net.address($listener)` to discover which port that was.
# Under `jennifer` (TinyGo build) every net call surfaces a
# friendly "not available" error - use `jennifer-go` for this.

use io;
use net;
use task;
use convert;

# ---- start the server ----
def listener as net.Listener init net.listen("127.0.0.1:0");
def addr as string init net.address($listener);

def server as task of string init spawn {
    def conn as net.Conn init net.accept($listener);
    def msg as bytes init net.readBytes($conn, 32);
    net.writeBytes($conn, $msg);
    net.close($conn);
    return convert.stringFromBytes($msg, "utf-8");
};

# ---- send + receive ----
def client as net.Conn init net.connect($addr);
net.writeBytes($client, convert.bytesFromString("hello, net", "utf-8"));
def reply as bytes init net.readBytes($client, 10);
net.close($client);

# ---- wait for the server side to finish so we can print its view ----
def seen as string init task.wait($server);
net.close($listener);

# ---- deterministic output (no port numbers) ----
io.printf("client received: %s\n", convert.stringFromBytes($reply, "utf-8"));
io.printf("server received: %s\n", $seen);
