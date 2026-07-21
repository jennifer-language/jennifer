#!/usr/bin/env -S jennifer run
# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 mplx <jennifer@mplx.dev>

/**
 * The amqp module (modules/amqp.j): an AMQP 0-9-1 client for RabbitMQ. Connect,
 * declare a queue, publish a few messages, then pull them back with Basic.Get
 * and acknowledge each. Needs the default `jennifer` binary (net) and a running
 * broker (default localhost:5672, guest/guest); pass host / user / password as
 * the first three arguments. Without a broker the connect throws, which this
 * demo catches and reports.
 * Run: jennifer run examples/modules/amqp_demo.j [host] [user] [password]
 * @module amqp_demo
 */
use io;
use os;
use convert;
import "../../modules/amqp.j" as amqp;

def host as string init "localhost";
def user as string init "guest";
def password as string init "guest";
if (len(os.ARGS) > 1) { $host = os.ARGS[1]; }
if (len(os.ARGS) > 2) { $user = os.ARGS[2]; }
if (len(os.ARGS) > 3) { $password = os.ARGS[3]; }

io.printf("connecting to %s (user %s) ...\n", $host, $user);
try {
    def c as amqp.Conn init amqp.connect(amqp.options($host, $user, $password));
    io.printf("connected; handshake ok\n");

    def qi as amqp.QueueInfo init amqp.declareQueue($c, "jennifer.demo", false);
    io.printf("queue %s: %d ready, %d consumers\n", $qi.name, $qi.messageCount, $qi.consumerCount);

    def payloads as list of string init ["one", "two", "three"];
    for (def p in $payloads) {
        amqp.publishText($c, "", "jennifer.demo", $p);
    }
    io.printf("published %d messages\n", len($payloads));

    def more as bool init true;
    def count as int init 0;
    repeat {
        def m as amqp.Message init amqp.get($c, "jennifer.demo", false);
        if ($m.empty) {
            $more = false;
        } else {
            io.printf("  got [tag %d] %s\n", $m.deliveryTag, convert.stringFromBytes($m.body, "utf-8"));
            amqp.ack($c, $m.deliveryTag);
            $count = $count + 1;
        }
    } until (not $more);
    io.printf("consumed %d messages\n", $count);

    amqp.close($c);
    io.printf("closed\n");
} catch (e) {
    io.printf("amqp unavailable: %s\n", $e.message);
}
