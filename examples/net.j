# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 <developer@mplx.eu>
#
# net.j - exercises the `net` library. A tiny in-process TCP echo: a
# `spawn`'d server accepts one connection and echoes whatever it reads;
# the main flow connects, sends a message, reads it back, and prints it.
# Then a short tour of the TLS client surface (connectTLS / startTLS /
# net.TLSOptions).
#
# Uses `:0` on the server so the kernel picks an ephemeral port, then
# `net.address($listener)` to discover which port that was. Under
# `jennifer-tiny` (TinyGo build) every net call surfaces a friendly
# "not available" error - use the default `jennifer` for this.

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

# ---- TLS client surface ----
# There is no in-process TLS server (net.listen is plaintext), so a real
# handshake talks to an external server. The client shape looks like:
#
#   def c as net.Conn init net.connectTLS("smtp.example.com:465");   # implicit TLS
#   net.writeBytes($c, ...); net.readBytes($c, n); net.close($c);    # same as plaintext
#
#   # STARTTLS: upgrade an open plaintext connection in place; the host
#   # is reused from connect(), so it isn't repeated:
#   def p as net.Conn init net.connect("smtp.example.com:587");
#   # ... EHLO / STARTTLS / read the "220 ready" line ...
#   $p = net.startTLS($p);
#
#   # net.TLSOptions (optional trailing arg): verification is on by
#   # default. Trust a self-signed cert with a PEM caCert (the secure
#   # path), or skipVerify to accept any cert (dev / testing only):
#   def o as net.TLSOptions;                # zero: skipVerify false, caCert empty
#   $o.caCert = fs.readBytes("server-ca.pem");
#   def s as net.Conn init net.connectTLS("localhost:8443", $o);
#
# The calls below just show the strict validation, which runs offline:

try {
    net.connectTLS("localhost");   # no port
    io.printf("missing port: not rejected\n");
} catch (portErr) {
    io.printf("connectTLS rejects a missing port\n");
}

def badca as net.TLSOptions;
$badca.caCert = convert.bytesFromString("not a certificate", "utf-8");
try {
    net.connectTLS("localhost:8443", $badca);
    io.printf("bad caCert: not rejected\n");
} catch (caErr) {
    io.printf("connectTLS rejects an invalid caCert PEM\n");
}
