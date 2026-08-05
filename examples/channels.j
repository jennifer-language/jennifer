# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

# CSP channels: streaming values between goroutines with `spawn` + `channel`.

use io;
use channel;
use task;

# 1. Producer / consumer over an unbuffered channel. The producer sends five
# values and closes; the consumer drains until the close raises, then stops.
def ch as channel of int init channel.make(0);
def producer as task of int init spawn {
    def i as int init 0;
    while ($i < 5) {
        channel.send($ch, $i * 10);
        $i = $i + 1;
    }
    channel.close($ch);
    return 0;
};
def sum as int init 0;
try {
    while (true) {
        $sum = $sum + channel.recv($ch);
    }
} catch (e) {
    # channel closed and drained
}
task.wait($producer);
io.printf("producer/consumer sum = %d\n", $sum);

# 2. Value semantics: the value is copied at send, so a later mutation by the
# sender does not reach the receiver.
def box as channel of list of int init channel.make(1);
def xs as list of int init [1, 2, 3];
channel.send($box, $xs);
$xs[0] = 999;
def got as list of int init channel.recv($box);
io.printf("sent copy = %v (sender mutated to %v)\n", $got, $xs);

# 3. Fan-in with select: merge two producers into one consumer.
def a as channel of int init channel.make(0);
def b as channel of int init channel.make(0);
def pa as task of int init spawn { channel.send($a, 100); channel.close($a); return 0; };
def pb as task of int init spawn { channel.send($b, 200); channel.close($b); return 0; };
def total as int init 0;
try {
    while (true) {
        $total = $total + channel.select([$a, $b]);
    }
} catch (e) {
    # both inputs closed
}
task.wait($pa);
task.wait($pb);
io.printf("fan-in select total = %d\n", $total);
