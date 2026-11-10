#!/usr/bin/env -S jennifer run
# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <mplx@jennifer-lang.dev>

/**
 * Request / reply and publish / subscribe with the zmq module, both ends in one
 * program over loopback: a spawned task serves, the main flow is the client.
 * The same sockets talk to libzmq, pyzmq or any other ZMTP 3 peer; point
 * `connect` at its "host:port". Needs the default `jennifer` binary (the module
 * uses `net`).
 * @module zmq_demo
 */
use io;
use convert;
use task;
use strings;
import "../../modules/zmq.j" as zmq;

func b(s as string) {
    return convert.bytesFromString("utf-8", $s);
}

# --- REQ / REP: a tiny upper-casing service ------------------------------------

def l as zmq.Listener init zmq.listen("REP", "127.0.0.1:0");
def addr as string init zmq.address($l);
def server as task of null init spawn {
    def s as zmq.Socket init zmq.accept($l);
    for (def i in 0..2) {
        def req as zmq.Message init zmq.receive($s);
        zmq.reply($s, $req, [b(strings.upper(zmq.text($req)))]);
    }
    zmq.close($s);
    return;
};

def c as zmq.Socket init zmq.connect("REQ", $addr);
for (def word in ["hello", "zeromq"]) {
    zmq.sendText($c, $word);
    io.printf("%s -> %s\n", $word, zmq.text(zmq.receive($c)));
}
zmq.close($c);
task.wait($server);
zmq.closeListener($l);

# --- PUB / SUB: topic frame + body frame ------------------------------------------

def pl as zmq.Listener init zmq.listen("PUB", "127.0.0.1:0");
def paddr as string init zmq.address($pl);
def subscriber as task of list of string init spawn {
    def s as zmq.Socket init zmq.subscribe(zmq.connect("SUB", $paddr), "sensor.");
    def got as list of string init [];
    for (def i in 0..2) {
        def m as zmq.Message init zmq.receive($s);
        $got[] = zmq.text($m) + " = " + convert.stringFromBytes("utf-8", $m.frames[1]);
    }
    zmq.close($s);
    return $got;
};
def pub as zmq.Socket init zmq.accept($pl);
zmq.send($pub, [b("log.debug"), b("not subscribed")]);
zmq.send($pub, [b("sensor.temp"), b("21.5")]);
zmq.send($pub, [b("sensor.humidity"), b("40")]);
for (def line in task.wait($subscriber)) {
    io.printf("%s\n", $line);
}
zmq.close($pub);
zmq.closeListener($pl);
